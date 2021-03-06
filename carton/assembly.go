/*
** Copyright [2013-2017] [Megam Systems]
**
** Licensed under the Apache License, Version 2.0 (the "License");
** you may not use this file except in compliance with the License.
** You may obtain a copy of the License at
**
** http://www.apache.org/licenses/LICENSE-2.0
**
** Unless required by applicable law or agreed to in writing, software
** distributed under the License is distributed on an "AS IS" BASIS,
** WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
** See the License for the specific language governing permissions and
** limitations under the License.
 */
package carton

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/virtengine/libgo/api"
	"github.com/virtengine/libgo/events"
	"github.com/virtengine/libgo/events/alerts"
	"github.com/virtengine/libgo/pairs"
	"github.com/virtengine/libgo/utils"
	constants "github.com/virtengine/libgo/utils"
	lb "github.com/virtengine/vertice/logbox"
	"github.com/virtengine/vertice/meta"
	"github.com/virtengine/vertice/provision"
	"gopkg.in/yaml.v2"
	// "github.com/virtengine/libgo/cmd"
)

const (
	ASSEMBLYBUCKET        = "assembly"
	ASM_UPDATE            = "/assembly/update"
	SSHKEY                = "sshkey"
	VNCPORT               = "vncport"
	VNCHOST               = "vnchost"
	INSTANCE_ID           = "instance_id"
	INSTANCE_PORTS        = "instance_ports"
	BACKUP                = "backup"
	YES                   = "yes"
	REGION                = "region"
	QUOTAID               = "quota_id"
	FLAVOR_ID             = "flavor_id"
	VM_CPU_COST           = "vm_cpu_cost_per_hour"
	VM_MEMORY_COST        = "vm_ram_cost_per_hour"
	VM_DISK_COST          = "vm_disk_cost_per_hour"
	CONTAINER_CPU_COST    = "container_cpu_cost_per_hour"
	CONTAINER_MEMORY_COST = "container_memory_cost_per_hour"
	CONTAINER_DISK_COST   = "container_disk_cost_per_hour"
)

type Policy struct {
	Name       string          `json:"name"`
	Type       string          `json:"ptype"`
	Resources  pairs.JsonPairs `json:"resources"`
	Rules      pairs.JsonPairs `json:"rules"`
	Properties pairs.JsonPairs `json:"properties"`
	Status     string          `json:"status"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

type Assembly struct {
	Id           string                `json:"id" cql:"id"`
	OrgId        string                `json:"org_id" cql:"org_id"`
	AccountId    string                `json:"account_id" cql:"account_id"`
	Name         string                `json:"name" cql:"name"`
	JsonClaz     string                `json:"json_claz" cql:"json_claz"`
	Tosca        string                `json:"tosca_type" cql:"tosca_type"`
	Status       string                `json:"status" cql:"status"`
	State        string                `json:"state" cql:"state"`
	CreatedAt    string                `json:"created_at" cql:"created_at"`
	Inputs       pairs.JsonPairs       `json:"inputs" cql:"inputs"`
	Outputs      pairs.JsonPairs       `json:"outputs" cql:"outputs"`
	Policies     []*Policy             `json:"policies" cql:"-"`
	ComponentIds []string              `json:"components" cql:"components"`
	Components   map[string]*Component `json:"-" cql:"-"`
}

type ApiAssembly struct {
	JsonClaz string     `json:"json_claz"`
	Results  []Assembly `json:"results"`
}

func (a *Assembly) String() string {
	if d, err := yaml.Marshal(a); err != nil {
		return err.Error()
	} else {
		return string(d)
	}
}

func get(args api.ApiArgs, ay string) (*Assembly, error) {
	cl := api.NewClient(args, "/assembly/"+ay)
	response, err := cl.Get()
	if err != nil {
		return nil, err
	}

	ac := &ApiAssembly{}
	//log.Debugf("Response %s :  (%s)",cmd.Colorfy("[Body]", "green", "", "bold"),string(htmlData))
	err = json.Unmarshal(response, ac)
	if err != nil {
		return nil, err
	}
	a := ac.Results[0]
	return a.dig()
}

// get all records in the assembly table with master credentials
func (a *Assembly) gets(args api.ApiArgs) ([]Assembly, error) {
	cl := api.NewClient(args, "/assembly")
	response, err := cl.Get()
	if err != nil {
		return nil, err
	}
	ac := &ApiAssembly{}
	err = json.Unmarshal(response, ac)
	if err != nil {
		return nil, err
	}
	if len(ac.Results) > 0 {
		return ac.Results, nil
	}
	return nil, fmt.Errorf("No records found")
}

func (a *Assembly) dig() (*Assembly, error) {
	a.Components = make(map[string]*Component)
	for _, cid := range a.ComponentIds {
		if len(strings.TrimSpace(cid)) > 1 {
			if comp, err := NewComponent(cid, a.AccountId, a.OrgId); err != nil {
				log.Errorf("Failed to get component %s from scylla: %s.", cid, err.Error())
				return a, err
			} else {
				a.Components[cid] = comp
			}
		}
	}
	return a, nil
}

func (a *Assembly) update() error {
	args := newArgs(a.AccountId, a.OrgId)
	cl := api.NewClient(args, "/assembly/update")
	_, err := cl.Post(a)
	if err != nil {
		return err
	}
	return nil
}

func NewArgs(email, org string) api.ApiArgs {
	return newArgs(email, org)
}

func newArgs(email, org string) api.ApiArgs {
	return api.ApiArgs{
		Master_Key: meta.MC.MasterKey,
		Url:        meta.MC.Api,
		Email:      email,
		Org_Id:     org,
	}
}

//Assembly into a carton.
//a carton comprises of self contained boxes
func mkCarton(aies, ay, email string) (*Carton, error) {
	args := newArgs(email, "")
	act, err := new(Account).get(args)
	if err != nil {
		return nil, err
	}

	a, err := get(args, ay)
	if err != nil {
		return nil, err
	}

	args.Api_Key = act.ApiKey
	args.Org_Id = a.OrgId
	b, err := a.mkBoxes(aies, args)
	if err != nil {
		return nil, err
	}

	c := &Carton{
		Id:           ay,   //assembly id
		CartonsId:    aies, //assemblies id
		OrgId:        a.OrgId,
		Name:         a.Name,
		Tosca:        a.Tosca,
		AccountId:    a.AccountId,
		Authority:    act.States.Authority,
		ApiArgs:      args,
		ImageVersion: a.imageVersion(),
		DomainName:   a.domain(),
		SSH:          a.newSSH(),
		Provider:     a.provider(),
		PublicIp:     a.publicIp(),
		Region:       a.region(),
		Vnets:        a.vnets(),
		InstanceId:   a.instanceId(),
		PolicyOps:    a.policyOps(),
		Backup:       a.isBackup(),
		ImageName:    a.imageName(),
		StorageType:  a.storageType(),
		QuotaId:      a.quotaID(),
		Boxes:        &b,
		Status:       utils.Status(a.Status),
		State:        utils.State(a.State),
	}
	if len(a.flavorId()) > 0 {
		comp, err := a.newCompute()
		if err != nil {
			return nil, err
		}
		c.Compute = comp
	} else {
		c.Compute = a.compute()
	}
	return c, nil
}

//lets make boxes with components to be mutated later or, and the required
//information for a launch.
//A "colored component" externalized with what we need.
func (a *Assembly) mkBoxes(aies string, args api.ApiArgs) ([]provision.Box, error) {
	vnet := a.vnets()
	instanceId := a.instanceId()
	newBoxs := make([]provision.Box, 0, len(a.Components))
	for _, comp := range a.Components {
		if len(strings.TrimSpace(comp.Id)) > 1 {
			if b, err := comp.mkBox(); err != nil {
				return nil, err
			} else {
				b.CartonId = a.Id
				b.CartonsId = aies
				b.CartonName = a.Name
				b.AccountId = a.AccountId
				b.OrgId = a.OrgId
				b.ApiArgs = args
				b.StorageType = a.storageType()
				if len(strings.TrimSpace(b.Provider)) <= 0 {
					b.Provider = a.provider()
				}
				if len(strings.TrimSpace(b.PublicIp)) <= 0 {
					b.PublicIp = a.publicIp()
				}
				if b.Repo.IsEnabled() {
					b.Repo.Hook.CartonId = a.Id //this is screwy, why do we need it.
					b.Repo.Hook.BoxId = comp.Id
				}
				if len(a.flavorId()) > 0 {
					c, err := a.newCompute()
					if err != nil {
						return nil, err
					}
					b.Compute = c
				} else {
					b.Compute = a.compute()
				}
				b.PolicyOps = a.policyOps()
				b.SSH = a.newSSH()
				b.Region = a.region()
				b.Status = utils.Status(a.Status)
				b.State = utils.State(a.State)
				b.Vnets = vnet
				b.InstanceId = instanceId
				b.QuotaId = a.quotaID()
				newBoxs = append(newBoxs, b)
			}
		}
	}
	return newBoxs, nil
}

//Temporary hack to create an assembly from its id.
//This is used by SetStatus.
//We need add a Notifier interface duck typed by Box and Carton ?
func NewAssembly(id, email, org string) (*Assembly, error) {
	args := newArgs(email, org)
	return get(args, id)
}

func AssemblyBox() ([]Assembly, error) {
	return new(Assembly).gets(newArgs(meta.MC.MasterUser, ""))
}

func NewCarton(aies, ay, email string) (*Carton, error) {
	return mkCarton(aies, ay, email)
}

func (a *Assembly) NukeKeysInputs(m string) error {
	if len(m) > 0 {
		log.Debugf("nuke keys from inputs in cassandra [%s]", m)
		a.Inputs.NukeKeys(m) //just nuke the matching output key:
		return a.update()
	}
	return provision.ErrNoOutputsFound
}

func (a *Assembly) SetStatus(status utils.Status) error {
	LastStatusUpdate := time.Now().Local().Format(time.RFC822)
	m := make(map[string][]string, 2)
	m["lastsuccessstatusupdate"] = []string{LastStatusUpdate}
	m["status"] = []string{status.String()}
	a.Inputs.NukeAndSet(m) //just nuke the matching output key:
	a.Status = status.String()
	err := a.update()
	if err != nil {
		return err
	}
	return a.trigger_event(status)
}

func (a *Assembly) SetState(state utils.State) error {
	a.State = state.String()
	return a.update()
}

func (a *Assembly) Trigger_event(status utils.Status) error {
	return a.trigger_event(status)
}

func (a *Assembly) trigger_event(status utils.Status) error {
	mi := make(map[string]string)
	js := make(pairs.JsonPairs, 0)
	m := make(map[string][]string, 2)
	m["status"] = []string{status.String()}
	m["description"] = []string{status.Description(a.Name)}
	js.NukeAndSet(m) //just nuke the matching output key:

	mi[constants.ASSEMBLY_ID] = a.Id
	mi[constants.ACCOUNT_ID] = a.AccountId
	mi[constants.EVENT_TYPE] = status.Event_type()

	newEvent := events.NewMulti(
		[]*events.Event{
			&events.Event{
				AccountsId:  a.AccountId,
				EventAction: alerts.STATUS,
				EventType:   constants.EventUser,
				EventData:   alerts.EventData{M: mi, D: js.ToString()},
				Timestamp:   time.Now().Local(),
			},
		})

	return newEvent.Write()
}

func DoneNotify(box *provision.Box, w io.Writer, evtAction alerts.EventAction, message string) error {
	fmt.Fprintf(w, lb.W(lb.DEPLOY, lb.INFO, fmt.Sprintf("--- done %s box ", box.GetFullName())))
	mi := make(map[string]string)
	mi[constants.VERTNAME] = box.GetFullName()
	mi[constants.VERTTYPE] = box.Tosca
	mi[constants.EMAIL] = box.AccountId
	if message != "" {
		mi[constants.ALERT_MESSAGE] = message
	}
	if evtAction == alerts.RUNNING {
		if box.SSH.Password != "" {
			pwd, _ := b64.StdEncoding.DecodeString(box.SSH.Password)
			mi[constants.INSTANCE_PASSWORD] = string(pwd)
		} else {
			mi[constants.SSH_KEY] = box.SSH.Prefix
		}
	}
	newEvent := events.NewMulti(
		[]*events.Event{
			&events.Event{
				AccountsId:  box.AccountId,
				EventAction: evtAction,
				EventType:   constants.EventMachine,
				EventData:   alerts.EventData{M: mi},
				Timestamp:   time.Now().Local(),
			},
		})
	fmt.Fprintf(w, lb.W(lb.DEPLOY, lb.INFO, fmt.Sprintf("--- done %s box OK", box.GetFullName())))
	return newEvent.Write()
}

//update outputs in scylla, nuke the matching keys available
func (a *Assembly) NukeAndSetOutputs(m map[string][]string) error {
	if len(m) > 0 {
		log.Debugf("nuke and set outputs in scylla [%s]", m)
		a.Outputs.NukeAndSet(m) //just nuke the matching output key:
		err := a.update()
		if err != nil {
			return err
		}
	} else {
		return provision.ErrNoOutputsFound
	}
	return nil
}

func (a *Assembly) Delete(asmid string) error {
	args := newArgs(a.AccountId, a.OrgId)
	cl := api.NewClient(args, "/assembly/"+asmid)
	_, err := cl.Delete()
	if err != nil {
		return err
	}
	return nil
}

func (a *Assembly) sshkey() string {
	return a.Inputs.Match(SSHKEY)
}

func (a *Assembly) domain() string {
	return a.Inputs.Match(DOMAIN)
}

func (a *Assembly) provider() string {
	return a.Inputs.Match(utils.PROVIDER)
}

func (a *Assembly) region() string {
	return a.Inputs.Match(REGION)
}

func (a *Assembly) vnets() map[string]string {
	v := make(map[string]string)
	v[utils.PUBLICIPV4] = a.ipv4Pub()
	v[utils.PRIVATEIPV4] = a.ipv4Pri()
	v[utils.PUBLICIPV6] = a.ipv6Pub()
	v[utils.PRIVATEIPV6] = a.ipv6Pri()
	return v
}

func (a *Assembly) ipv4Pub() string {
	return a.Inputs.Match(utils.PUBLICIPV4)
}

func (a *Assembly) ipv4Pri() string {
	return a.Inputs.Match(utils.PRIVATEIPV4)
}

func (a *Assembly) ipv6Pri() string {
	return a.Inputs.Match(utils.PRIVATEIPV6)
}

func (a *Assembly) ipv6Pub() string {
	return a.Inputs.Match(utils.PUBLICIPV6)
}

func (a *Assembly) publicIp() string {
	return a.Outputs.Match(utils.PUBLICIPV4)
}
func (a *Assembly) vncHost() string {
	return a.Outputs.Match(VNCHOST)
}
func (a *Assembly) vncPort() string {
	return a.Outputs.Match(VNCPORT)
}
func (a *Assembly) instanceId() string {
	return a.Outputs.Match(INSTANCE_ID)
}
func (a *Assembly) imageVersion() string {
	return a.Inputs.Match(IMAGE_VERSION)
}

func (a *Assembly) imageName() string {
	return a.Inputs.Match(BACKUPNAME)
}

func (a *Assembly) quotaID() string {
	return strings.TrimSpace(a.Inputs.Match(QUOTAID))
}

func (a *Assembly) QuotaId() string {
	return a.quotaID()
}

func (a *Assembly) IsQuota() bool {
	return len(a.quotaID()) > 0
}

func (a *Assembly) storageType() string {
	return strings.ToLower(a.Inputs.Match(utils.STORAGE_TYPE))
}

func (a *Assembly) isBackup() bool {
	return (strings.TrimSpace(a.Inputs.Match(BACKUP)) == YES)
}

func (a *Assembly) flavorId() string {
	return strings.TrimSpace(a.Inputs.Match(FLAVOR_ID))
}

func (a *Assembly) FlavorId() string {
	return strings.TrimSpace(a.Inputs.Match(FLAVOR_ID))
}

func (a *Assembly) newCompute() (provision.BoxCompute, error) {
	comp := provision.BoxCompute{}
	f, err := GetFlavor(a.AccountId, a.flavorId())
	if err != nil {
		return comp, err
	}
	return f.compute(), nil
}

func (a *Assembly) compute() provision.BoxCompute {
	return provision.BoxCompute{
		Cpushare: a.getCpushare(),
		Memory:   a.getMemory(),
		Swap:     a.getSwap(),
		HDD:      a.getHDD(),
	}
}

func (a *Assembly) newSSH() provision.BoxSSH {
	return provision.BoxSSH{
		User:     meta.MC.User,
		Prefix:   a.sshkey(),
		Password: a.password(),
	}
}

func (a *Assembly) password() string {
	return a.Inputs.Match(constants.ROOT_PASSWORD)
}

func (a *Assembly) getCpushare() string {
	return a.Inputs.Match(provision.CPU)
}

func (a *Assembly) getMemory() string {
	return a.Inputs.Match(provision.RAM)
}

func (a *Assembly) getSwap() string {
	return ""
}

//The default HDD is 10. we should configure it in the vertice.conf
func (a *Assembly) getHDD() string {
	if len(strings.TrimSpace(a.Inputs.Match(provision.HDD))) <= 0 {
		return "10"
	}
	return a.Inputs.Match(provision.HDD)
}

func (a *Assembly) GetVMCpuCost() string {
	return a.Inputs.Match(VM_CPU_COST)
}

func (a *Assembly) GetVMMemoryCost() string {
	return a.Inputs.Match(VM_MEMORY_COST)
}

func (a *Assembly) GetVMHDDCost() string {
	return a.Inputs.Match(VM_DISK_COST)
}

func (a *Assembly) GetContainerCpuCost() string {
	return a.Inputs.Match(CONTAINER_CPU_COST)
}

func (a *Assembly) GetContainerMemoryCost() string {
	return a.Inputs.Match(CONTAINER_MEMORY_COST)
}

func (a *Assembly) HostName() string {
	return a.Outputs.Match(VNCHOST)
}

func (a *Assembly) GetFullName() string {
	domain := a.domain()
	if len(strings.TrimSpace(domain)) > 0 {
		return strings.Join([]string{a.Name, domain}, ".")
	}
	return a.Name
}

func parseStringToStruct(str string, data interface{}) error {
	if err := json.Unmarshal([]byte(str), data); err != nil {
		return err
	}
	return nil
}

func (a *Assembly) UpdatePolicyStatus(index int, status utils.Status) error {
	a.Policies[index].Status = status.String()
	return a.update()
}

func (a *Assembly) policyOps() *provision.PolicyOps {
	for i, policy := range a.Policies {
		if policy.Status == "initializing" {
			return &provision.PolicyOps{
				Type:       policy.Type,
				Operation:  policy.Name,
				Index:      i,
				Rules:      policy.rules(),
				Properties: policy.properties(),
			}
		}
	}
	return nil
}

func (p *Policy) rules() map[string]string {
	return p.Rules.ToMap()
}

func (p *Policy) properties() map[string]string {
	return p.Properties.ToMap()
}

func (a *Assembly) SetStatusErr(status utils.Status, causeof error) error {
	LastStatusUpdate := time.Now().Local().Format(time.RFC822)
	m := make(map[string][]string, 2)
	m["lastsuccessstatusupdate"] = []string{LastStatusUpdate}
	m["status"] = []string{status.String()}
	a.Inputs.NukeAndSet(m) //just nuke the matching output key:
	a.Status = status.String()
	err := a.update()
	if err != nil {
		return err
	}
	return a.trigger_error_event(status, causeof)
}

func (a *Assembly) trigger_error_event(status utils.Status, causeof error) error {
	mi := make(map[string]string)
	js := make(pairs.JsonPairs, 0)
	m := make(map[string][]string, 2)
	m["status"] = []string{status.String()}
	m["description"] = []string{status.Description(causeof.Error())}
	js.NukeAndSet(m) //just nuke the matching output key:

	mi[constants.ASSEMBLY_ID] = a.Id
	mi[constants.ACCOUNT_ID] = a.AccountId
	mi[constants.EVENT_TYPE] = status.Event_type()

	newEvent := events.NewMulti(
		[]*events.Event{
			&events.Event{
				AccountsId:  a.AccountId,
				EventAction: alerts.STATUS,
				EventType:   constants.EventUser,
				EventData:   alerts.EventData{M: mi, D: js.ToString()},
				Timestamp:   time.Now().Local(),
			},
		})

	return newEvent.Write()
}
func (a *Assembly) IsAlive() bool {
	return !a.isPending() && !a.isDestroyed()
}

func (a *Assembly) isPending() bool {
	return a.State == constants.PREDEPLOY_ERROR || a.State == constants.PARKED || a.State == constants.INITIALIZING
}

func (a *Assembly) isDestroyed() bool {
	return a.State == constants.DESTROYING || a.State == constants.DESTROYED
}

func (a *Assembly) isStopped() bool {
	return a.State == constants.STOPPED || a.State == constants.STOPPING
}

func (a *Assembly) isSuspended() bool {
	return a.State == constants.SUSPENDED || a.State == constants.SUSPENDING
}

func (a *Assembly) IsContainer() bool {
	return strings.Split(a.Tosca, ".")[1] == constants.CONTAINER
}

func (a *Assembly) IsTopedo() bool {
	return strings.Split(a.Tosca, ".")[1] == constants.TORPEDO
}
func (a *Assembly) Resources(flv *Flavor) map[string]string {
	box := &provision.Box{}
	if flv != nil {
		box.Compute = flv.compute()
		return a.billaleResource(map[string]string{
			constants.CPU:         strconv.FormatInt(int64(box.GetCpushare()), 10),
			constants.RAM:         strconv.FormatInt(int64(box.GetMemory()), 10),
			constants.STORAGE:     strconv.FormatInt(int64(box.GetHDD()), 10),
			constants.CPU_COST:    flv.GetCpuCost(),
			constants.MEMORY_COST: flv.GetMemoryCost(),
			constants.DISK_COST:   flv.GetHDDCost(),
		})
	}
	return a.resources()
}

func (a *Assembly) resources() map[string]string {
	box := &provision.Box{Compute: a.compute()}
	r := map[string]string{
		constants.CPU:     strconv.FormatInt(int64(box.GetCpushare()), 10),
		constants.RAM:     strconv.FormatInt(int64(box.GetMemory()), 10),
		constants.STORAGE: strconv.FormatInt(int64(box.GetHDD()), 10),
	}
	if a.IsContainer() {
		r[constants.CPU_COST] = a.GetContainerCpuCost()
		r[constants.MEMORY_COST] = a.GetContainerMemoryCost()
	} else {
		r[constants.CPU_COST] = a.GetVMCpuCost()
		r[constants.MEMORY_COST] = a.GetVMMemoryCost()
		r[constants.DISK_COST] = a.GetVMHDDCost()
	}
	return a.billaleResource(r)
}

func (a *Assembly) billaleResource(r map[string]string) map[string]string {
	if a.isStopped() || a.isSuspended() {
		r[constants.CPU_COST] = "0"
		r[constants.MEMORY_COST] = "0"
		r[constants.RESOURCES] = "storage"
	} else {
		r[constants.RESOURCES] = "cpu.ram.storage"
	}
	return r
}
