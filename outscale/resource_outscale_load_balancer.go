package outscale

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	oscgo "github.com/outscale/osc-sdk-go/v2"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/terraform-providers/terraform-provider-outscale/utils"
)

func lb_sg_schema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Computed: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"security_group_name": {
					Type:     schema.TypeString,
					Computed: true,
				},
				"security_group_account_id": {
					Type:     schema.TypeString,
					Computed: true,
				},
			},
		},
	}
}

func resourceOutscaleOAPILoadBalancer() *schema.Resource {
	return &schema.Resource{
		Create: resourceOutscaleOAPILoadBalancerCreate,
		Read:   resourceOutscaleOAPILoadBalancerRead,
		Update: resourceOutscaleOAPILoadBalancerUpdate,
		Delete: resourceOutscaleOAPILoadBalancerDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"subregion_names": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				ForceNew: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"load_balancer_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"load_balancer_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"security_groups": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"subnets": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"tags": tagsListOAPISchema2(false),

			"dns_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"access_log": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"is_enabled": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"osu_bucket_name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"osu_bucket_prefix": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"publication_interval": {
							Type:     schema.TypeInt,
							Computed: true,
						},
					},
				},
			},
			"health_check": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"healthy_threshold": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"unhealthy_threshold": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"path": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"check_interval": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"port": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"protocol": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"timeout": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"backend_vm_ids": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"listeners": {
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: lb_listener_schema(false),
				},
			},
			"source_security_group": lb_sg_schema(),
			"net_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"application_sticky_cookie_policies": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cookie_name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"policy_name": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"load_balancer_sticky_cookie_policies": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"policy_name": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"request_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func expandStringList(ifs []interface{}) *[]string {
	r := make([]string, len(ifs))

	for k, v := range ifs {
		r[k] = v.(string)
	}
	return &r
}

func expandSetStringList(ifs *schema.Set) *[]string {
	r := make([]string, ifs.Len())

	for k, v := range ifs.List() {
		r[k] = v.(string)
	}
	return &r
}

// Flattens an array of Listeners into a []map[string]interface{}
func flattenOAPIListeners(list *[]oscgo.Listener) []map[string]interface{} {
	if list == nil {
		return make([]map[string]interface{}, 0)
	}

	result := make([]map[string]interface{}, 0, len(*list))

	for _, i := range *list {
		listener := map[string]interface{}{
			"backend_port":           int(*i.BackendPort),
			"backend_protocol":       *i.BackendProtocol,
			"load_balancer_port":     int(*i.LoadBalancerPort),
			"load_balancer_protocol": *i.LoadBalancerProtocol,
		}
		if i.ServerCertificateId != nil {
			listener["server_certificate_id"] =
				*i.ServerCertificateId
		}
		listener["policy_names"] = flattenStringList(i.PolicyNames)
		result = append(result, listener)
	}
	return result
}

func expandListeners(configured []interface{}) ([]*oscgo.Listener, error) {
	listeners := make([]*oscgo.Listener, 0, len(configured))

	for _, lRaw := range configured {
		data := lRaw.(map[string]interface{})

		ip := int32(data["backend_port"].(int))
		lp := int32(data["load_balancer_port"].(int))
		bproto := data["backend_protocol"].(string)
		lproto := data["load_balancer_protocol"].(string)
		l := &oscgo.Listener{
			BackendPort:          &ip,
			BackendProtocol:      &bproto,
			LoadBalancerPort:     &lp,
			LoadBalancerProtocol: &lproto,
		}

		if v, ok := data["server_certificate_id"]; ok && v != "" {
			vs := v.(string)
			l.ServerCertificateId = &vs
		}

		var valid bool
		if l.ServerCertificateId != nil && *l.ServerCertificateId != "" {
			// validate the protocol is correct
			for _, p := range []string{"https", "ssl"} {
				if (strings.ToLower(*l.BackendProtocol) == p) ||
					(strings.ToLower(*l.LoadBalancerProtocol) == p) {
					valid = true
				}
			}
		} else {
			valid = true
		}

		if valid {
			listeners = append(listeners, l)
		} else {
			return nil, fmt.Errorf("[ERR] ELB Listener: server_certificate_id may be set only when protocol is 'https' or 'ssl'")
		}
	}

	return listeners, nil
}

func expandListenerForCreation(configured []interface{}) ([]oscgo.ListenerForCreation, error) {
	listeners := make([]oscgo.ListenerForCreation, 0, len(configured))

	for _, lRaw := range configured {
		data := lRaw.(map[string]interface{})

		ip := int32(data["backend_port"].(int))
		lp := int32(data["load_balancer_port"].(int))
		bproto := data["backend_protocol"].(string)
		lproto := data["load_balancer_protocol"].(string)
		l := oscgo.ListenerForCreation{
			BackendPort:          ip,
			BackendProtocol:      &bproto,
			LoadBalancerPort:     lp,
			LoadBalancerProtocol: lproto,
		}

		if v, ok := data["server_certificate_id"]; ok && v != "" {
			vs := v.(string)
			l.ServerCertificateId = &vs
		}

		var valid bool
		if l.ServerCertificateId != nil && *l.ServerCertificateId != "" {
			// validate the protocol is correct
			for _, p := range []string{"https", "ssl"} {
				if (strings.ToLower(*l.BackendProtocol) == p) ||
					(strings.ToLower(l.LoadBalancerProtocol) == p) {
					valid = true
				}
			}
		} else {
			valid = true
		}

		if valid {
			listeners = append(listeners, l)
		} else {
			return nil, fmt.Errorf("[ERR] ELB Listener: server_certificate_id may be set only when protocol is 'https' or 'ssl'")
		}
	}

	return listeners, nil
}

func mk_elem(computed bool, required bool, optional bool,
	t schema.ValueType) *schema.Schema {
	if computed {
		return &schema.Schema{
			Type:     t,
			Computed: true,
		}
	} else if required {
		return &schema.Schema{
			Type:     t,
			Required: true,
		}
	} else {
		return &schema.Schema{
			Type:     t,
			Optional: true,
		}
	}
}

func lb_listener_schema(computed bool) map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"backend_port": mk_elem(computed, !computed, false,
			schema.TypeInt),
		"backend_protocol": mk_elem(computed, !computed, false,
			schema.TypeString),
		"load_balancer_port": mk_elem(computed, !computed, false,
			schema.TypeInt),
		"load_balancer_protocol": mk_elem(computed, !computed, false,
			schema.TypeString),
		"server_certificate_id": mk_elem(computed, false, !computed,
			schema.TypeString),
		"policy_names": {
			Type:     schema.TypeList,
			Computed: true,
			Elem:     &schema.Schema{Type: schema.TypeString},
		},
	}
}

func resourceOutscaleOAPILoadBalancerCreate(d *schema.ResourceData, meta interface{}) error {
	return resourceOutscaleOAPILoadBalancerCreate_(d, meta, false)
}

func resourceOutscaleOAPILoadBalancerCreate_(d *schema.ResourceData, meta interface{}, isUpdate bool) error {
	conn := meta.(*OutscaleClient).OSCAPI

	req := &oscgo.CreateLoadBalancerRequest{}

	listeners, err := expandListenerForCreation(d.Get("listeners").(*schema.Set).List())
	if err != nil {
		return err
	}

	req.Listeners = listeners

	if v, ok := d.GetOk("load_balancer_name"); ok {
		req.LoadBalancerName = v.(string)
	}

	if v, ok := d.GetOk("tags"); ok {
		r := tagsFromSliceMap(v.(*schema.Set))
		req.Tags = &r
	}

	if v, ok := d.GetOk("load_balancer_type"); ok {
		s := v.(string)
		req.LoadBalancerType = &s
	}

	if v, ok := d.GetOk("security_groups"); ok {
		req.SecurityGroups = expandSetStringList(v.(*schema.Set))
	}

	v_sb, sb_ok := d.GetOk("subnets")
	if sb_ok {
		req.Subnets = expandStringList(v_sb.([]interface{}))
	}

	v_srn, srn_ok := d.GetOk("subregion_names")
	if sb_ok && srn_ok {
		return fmt.Errorf("can't use both 'subregion_names' and 'subnets'")
	}

	if srn_ok && sb_ok == false {
		req.SubregionNames = expandStringList(v_srn.([]interface{}))
	}

	log.Printf("[DEBUG] Load Balancer request configuration: %#v", *req)
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, _, err = conn.LoadBalancerApi.CreateLoadBalancer(
			context.Background()).
			CreateLoadBalancerRequest(*req).Execute()

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "CertificateNotFound") {
				return resource.RetryableError(
					fmt.Errorf("[WARN] Error creating Load Balancer Listener with SSL Cert, retrying: %s", err))
			}
			if strings.Contains(fmt.Sprint(err), "Throttling") {
				return resource.RetryableError(
					fmt.Errorf("[WARN] Error creating Load Balancer Listener with SSL Cert, retrying: %s", err))
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Assign the lbu's unique identifier for use later
	d.SetId(req.LoadBalancerName)
	log.Printf("[INFO] Load Balancer ID: %s", d.Id())

	if err := d.Set("listeners", make([]map[string]interface{}, 0)); err != nil {
		return err
	}

	return resourceOutscaleOAPILoadBalancerRead(d, meta)
}

func flattenStringList(list *[]string) []interface{} {
	if list == nil {
		return make([]interface{}, 0)
	}
	vs := make([]interface{}, 0, len(*list))
	for _, v := range *list {
		vs = append(vs, v)
	}
	return vs
}

func readResourceLb(conn *oscgo.APIClient, elbName string) (*oscgo.LoadBalancer, *oscgo.ReadLoadBalancersResponse, error) {
	filter := &oscgo.FiltersLoadBalancer{
		LoadBalancerNames: &[]string{elbName},
	}

	req := oscgo.ReadLoadBalancersRequest{
		Filters: filter,
	}

	var resp oscgo.ReadLoadBalancersResponse
	var err error
	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, _, err = conn.LoadBalancerApi.ReadLoadBalancers(
			context.Background()).
			ReadLoadBalancersRequest(req).Execute()
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "Throttling:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return nil, nil, fmt.Errorf("Error retrieving Load Balancer: %s", err)
	}

	if err := utils.IsResponseEmptyOrMutiple(len(resp.GetLoadBalancers()), "LoadBalancer"); err != nil {
		return nil, nil, err
	}

	lb := (*resp.LoadBalancers)[0]
	return &lb, &resp, nil
}

func resourceOutscaleOAPILoadBalancerRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI
	elbName := d.Id()

	lb, _, err := readResourceLb(conn, elbName)
	if err != nil {
		return err
	}

	d.Set("subregion_names", flattenStringList(lb.SubregionNames))
	d.Set("dns_name", lb.DnsName)
	d.Set("health_check", flattenOAPIHealthCheck(lb.HealthCheck))
	d.Set("access_log", flattenOAPIAccessLog(lb.AccessLog))

	d.Set("backend_vm_ids", flattenStringList(lb.BackendVmIds))
	if err := d.Set("listeners", flattenOAPIListeners(lb.Listeners)); err != nil {
		log.Printf("[DEBUG] out err %v", err)
		return err
	}
	d.Set("load_balancer_name", lb.LoadBalancerName)

	if lb.Tags != nil {
		ta := make([]map[string]interface{}, len(*lb.Tags))
		for k1, v1 := range *lb.Tags {
			t := make(map[string]interface{})
			t["key"] = v1.Key
			t["value"] = v1.Value
			ta[k1] = t
		}

		d.Set("tags", ta)
	} else {
		d.Set("tags", make([]map[string]interface{}, 0))

	}

	if lb.ApplicationStickyCookiePolicies != nil {
		app := make([]map[string]interface{},
			len(*lb.ApplicationStickyCookiePolicies))
		for k, v := range *lb.ApplicationStickyCookiePolicies {
			a := make(map[string]interface{})
			a["cookie_name"] = v.CookieName
			a["policy_name"] = v.PolicyName
			app[k] = a
		}
		d.Set("application_sticky_cookie_policies", app)
	}
	if lb.LoadBalancerStickyCookiePolicies != nil {
		lbc := make([]map[string]interface{},
			len(*lb.LoadBalancerStickyCookiePolicies))
		for k, v := range *lb.LoadBalancerStickyCookiePolicies {
			a := make(map[string]interface{})
			a["policy_name"] = v.PolicyName
			lbc[k] = a
		}
		d.Set("load_balancer_sticky_cookie_policies", lbc)
	}

	d.Set("load_balancer_type", lb.LoadBalancerType)
	if lb.SecurityGroups != nil {
		d.Set("security_groups", flattenStringList(lb.SecurityGroups))
	} else {
		d.Set("security_groups", make([]map[string]interface{}, 0))
	}
	ssg := make(map[string]string)
	if lb.SourceSecurityGroup != nil {
		ssg["security_group_name"] = *lb.SourceSecurityGroup.SecurityGroupName
		ssg["security_group_account_id"] = *lb.SourceSecurityGroup.SecurityGroupAccountId
	}
	d.Set("source_security_group", ssg)
	d.Set("subnets", flattenStringList(lb.Subnets))
	d.Set("net_id", lb.NetId)

	return nil
}

func resourceOutscaleOAPILoadBalancerUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI
	var err error
	d.Partial(true)

	if d.HasChange("security_groups") {
		req := oscgo.UpdateLoadBalancerRequest{
			LoadBalancerName: d.Id(),
		}
		nSg, _ := d.GetOk("security_groups")
		req.SecurityGroups = expandSetStringList(nSg.(*schema.Set))

		var err error
		err = resource.Retry(1*time.Minute, func() *resource.RetryError {
			_, _, err = conn.LoadBalancerApi.UpdateLoadBalancer(
				context.Background()).UpdateLoadBalancerRequest(req).Execute()
			if err != nil {
				if strings.Contains(err.Error(), "Throttling:") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("Failure updating SecurityGroups: %s", err)
		}
		d.SetPartial("security_groups")
	}

	if d.HasChange("tags") {
		oraw, nraw := d.GetChange("tags")
		o := oraw.(*schema.Set)
		n := nraw.(*schema.Set)
		create := tagsFromSliceMap(n)
		var remove []oscgo.ResourceLoadBalancerTag
		for _, t := range o.List() {
			tag := t.(map[string]interface{})
			s := tag["key"].(string)
			remove = append(remove,
				oscgo.ResourceLoadBalancerTag{
					Key: &s,
				})
		}
		if len(remove) < 1 {
			goto skip_delete
		}

		err = resource.Retry(60*time.Second, func() *resource.RetryError {
			_, _, err := conn.LoadBalancerApi.DeleteLoadBalancerTags(
				context.Background()).
				DeleteLoadBalancerTagsRequest(
					oscgo.DeleteLoadBalancerTagsRequest{
						LoadBalancerNames: []string{d.Id()},
						Tags:              remove,
					}).Execute()
			if err != nil {
				if strings.Contains(fmt.Sprint(err), ".NotFound") {
					return resource.RetryableError(err) // retry
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})
		if err != nil {
			return err
		}

	skip_delete:
		if len(create) < 1 {
			goto skip_create
		}

		err = resource.Retry(60*time.Second, func() *resource.RetryError {
			_, _, err := conn.LoadBalancerApi.CreateLoadBalancerTags(
				context.Background()).
				CreateLoadBalancerTagsRequest(
					oscgo.CreateLoadBalancerTagsRequest{
						LoadBalancerNames: []string{d.Id()},
						Tags:              create,
					}).Execute()
			if err != nil {
				if strings.Contains(fmt.Sprint(err), ".NotFound") {
					return resource.RetryableError(err) // retry
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})
		if err != nil {
			return err
		}

	skip_create:
	}

	if d.HasChange("listeners") {
		o, n := d.GetChange("listeners")
		os := o.(*schema.Set).List()
		ns := n.(*schema.Set).List()

		log.Printf("[DEBUG] it change !: %v %v", os, ns)
		remove, _ := expandListeners(os)
		add, _ := expandListenerForCreation(ns)

		if len(remove) > 0 {
			ports := make([]int32, 0, len(remove))
			for _, listener := range remove {
				ports = append(ports, *listener.LoadBalancerPort)
			}

			req := oscgo.DeleteLoadBalancerListenersRequest{
				LoadBalancerName:  d.Id(),
				LoadBalancerPorts: ports,
			}

			log.Printf("[DEBUG] Load Balancer Delete Listeners")

			var err error
			err = resource.Retry(5*time.Minute, func() *resource.RetryError {
				_, _, err = conn.ListenerApi.DeleteLoadBalancerListeners(
					context.Background()).
					DeleteLoadBalancerListenersRequest(req).
					Execute()

				if err != nil {
					if strings.Contains(err.Error(), "Throttling:") {
						return resource.RetryableError(err)
					}
					return resource.NonRetryableError(err)
				}
				return nil
			})

			if err != nil {
				return fmt.Errorf("Failure removing outdated Load Balancer listeners: %s", err)
			}
		}

		if len(add) > 0 {
			req := oscgo.CreateLoadBalancerListenersRequest{
				LoadBalancerName: d.Id(),
				Listeners:        add,
			}

			// Occasionally AWS will error with a 'duplicate listener', without any
			// other listeners on the Load Balancer. Retry here to eliminate that.
			var err error
			err = resource.Retry(5*time.Minute, func() *resource.RetryError {
				log.Printf("[DEBUG] Load Balancer Create Listeners")
				_, _, err = conn.ListenerApi.CreateLoadBalancerListeners(
					context.Background()).CreateLoadBalancerListenersRequest(req).Execute()
				if err != nil {
					if strings.Contains(fmt.Sprint(err), "DuplicateListener") {
						log.Printf("[DEBUG] Duplicate listener found for ELB (%s), retrying", d.Id())
						return resource.RetryableError(err)
					}
					if strings.Contains(fmt.Sprint(err), "CertificateNotFound") && strings.Contains(fmt.Sprint(err), "Server Certificate not found for the key: arn") {
						log.Printf("[DEBUG] SSL Cert not found for given ARN, retrying")
						return resource.RetryableError(err)
					}
					if strings.Contains(fmt.Sprint(err), "Throttling") {
						return resource.RetryableError(
							fmt.Errorf("[WARN] Error creating ELB Listener with SSL Cert, retrying: %s", err))
					}
					return resource.NonRetryableError(err)
				}
				// Successful creation
				return nil
			})
			if err != nil {
				return fmt.Errorf("Failure adding new or updated Load Balancer listeners: %s", err)
			}
		}

		d.SetPartial("listeners")
	}

	if d.HasChange("backend_vm_ids") {
		o, n := d.GetChange("backend_vm_ids")
		os := o.(*schema.Set)
		ns := n.(*schema.Set)
		remove := expandInstanceString(os.Difference(ns).List())
		add := expandInstanceString(ns.Difference(os).List())

		if len(add) > 0 {

			req := oscgo.RegisterVmsInLoadBalancerRequest{
				LoadBalancerName: d.Id(),
				BackendVmIds:     add,
			}

			var err error
			err = resource.Retry(5*time.Minute, func() *resource.RetryError {
				_, _, err = conn.LoadBalancerApi.
					RegisterVmsInLoadBalancer(context.Background()).
					RegisterVmsInLoadBalancerRequest(req).
					Execute()

				if err != nil {
					if strings.Contains(fmt.Sprint(err), "Throttling") {
						return resource.RetryableError(
							fmt.Errorf("[WARN] Error, retrying: %s", err))
					}
					return resource.NonRetryableError(err)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("Failure registering instances with Load Balancer: %s", err)
			}
		}
		if len(remove) > 0 {
			req := oscgo.DeregisterVmsInLoadBalancerRequest{
				LoadBalancerName: d.Id(),
				BackendVmIds:     remove,
			}

			var err error
			err = resource.Retry(5*time.Minute, func() *resource.RetryError {
				_, _, err := conn.LoadBalancerApi.
					DeregisterVmsInLoadBalancer(
						context.Background()).
					DeregisterVmsInLoadBalancerRequest(req).
					Execute()

				if err != nil {
					if strings.Contains(err.Error(), "Throttling:") {
						return resource.RetryableError(err)
					}
					return resource.NonRetryableError(err)
				}
				return nil
			})

			if err != nil {
				return fmt.Errorf("Failure deregistering instances from Load Balancer: %s", err)
			}
		}

		d.SetPartial("backend_vm_ids")
	}

	if d.HasChange("health_check") {
		hc := d.Get("health_check").([]interface{})
		if len(hc) > 0 {
			check := hc[0].(map[string]interface{})
			req := oscgo.UpdateLoadBalancerRequest{
				LoadBalancerName: d.Id(),
				HealthCheck: &oscgo.HealthCheck{
					HealthyThreshold:   int32(check["healthy_threshold"].(int)),
					UnhealthyThreshold: int32(check["unhealthy_threshold"].(int)),
					CheckInterval:      int32(check["check_interval"].(int)),
					Protocol:           check["protocol"].(string),
					Port:               int32(check["port"].(int)),
					Timeout:            int32(check["timeout"].(int)),
				},
			}
			if check["path"] != nil {
				p := check["path"].(string)
				req.HealthCheck.Path = &p
			}

			var err error

			err = resource.Retry(5*time.Minute, func() *resource.RetryError {
				_, _, err = conn.LoadBalancerApi.UpdateLoadBalancer(
					context.Background()).UpdateLoadBalancerRequest(req).
					Execute()

				if err != nil {
					if strings.Contains(err.Error(), "Throttling:") {
						return resource.RetryableError(err)
					}
					return resource.NonRetryableError(err)
				}
				return nil
			})

			if err != nil {
				return fmt.Errorf("Failure configuring health check for Load Balancer: %s", err)
			}
			d.SetPartial("health_check")
		}
	}

	if d.HasChange("access_log") {
		acg := d.Get("access_log").([]interface{})
		if len(acg) > 0 {

			aclg := acg[0].(map[string]interface{})
			isEnabled := aclg["is_enabled"].(bool)
			osuBucketName := aclg["osu_bucket_name"].(string)
			osuBucketPrefix := aclg["osu_bucket_prefix"].(string)
			publicationInterval := int32(aclg["publication_interval"].(int))
			req := oscgo.UpdateLoadBalancerRequest{
				LoadBalancerName: d.Id(),
				AccessLog: &oscgo.AccessLog{
					IsEnabled:           &isEnabled,
					OsuBucketName:       &osuBucketName,
					OsuBucketPrefix:     &osuBucketPrefix,
					PublicationInterval: &publicationInterval,
				},
			}

			var err error

			err = resource.Retry(5*time.Minute, func() *resource.RetryError {
				_, _, err = conn.LoadBalancerApi.UpdateLoadBalancer(
					context.Background()).UpdateLoadBalancerRequest(req).Execute()

				if err != nil {
					if strings.Contains(err.Error(), "Throttling:") {
						return resource.RetryableError(err)
					}
					return resource.NonRetryableError(err)
				}
				return nil
			})

			if err != nil {
				return fmt.Errorf("Failure configuring access log for Load Balancer: %s", err)
			}
			d.SetPartial("access_log")
		}
	}

	d.SetPartial("listeners")
	d.SetPartial("application_sticky_cookie_policies")
	d.SetPartial("load_balancer_sticky_cookie_policies")

	d.Partial(false)

	return resourceOutscaleOAPILoadBalancerRead(d, meta)
}

func resourceOutscaleOAPILoadBalancerDelete(d *schema.ResourceData, meta interface{}) error {
	return resourceOutscaleOAPILoadBalancerDelete_(d, meta, true)
}

func resourceOutscaleOAPILoadBalancerDelete_(d *schema.ResourceData, meta interface{}, needupdate bool) error {
	conn := meta.(*OutscaleClient).OSCAPI

	log.Printf("[INFO] Deleting Load Balancer: %s", d.Id())

	// Destroy the load balancer
	req := oscgo.DeleteLoadBalancerRequest{
		LoadBalancerName: d.Id(),
	}

	var err error

	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, _, err = conn.LoadBalancerApi.DeleteLoadBalancer(
			context.Background()).DeleteLoadBalancerRequest(req).Execute()
		if err != nil {
			if strings.Contains(err.Error(), "Throttling:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error deleting Load Balancer: %s", err)
	}

	if needupdate {
		d.SetId("")
	}

	stateConf := &resource.StateChangeConf{
		Pending: []string{"ready"},
		Target:  []string{},
		Refresh: func() (interface{}, string, error) {
			lb, _, _ := readResourceLb(conn, d.Id())
			if lb == nil {
				return nil, "", nil
			}
			return lb, "ready", nil
		},
		Timeout: 5 * time.Minute,
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf("Error waiting for load balancer (%s) to become null: %s", d.Id(), err)
	}

	return nil
}

// Expands an array of String Instance IDs into a []Instances
func expandInstanceString(list []interface{}) []string {
	result := make([]string, 0, len(list))
	for _, i := range list {
		result = append(result, i.(string))
	}
	return result
}

func formatInt32(n int32) string {
	return strconv.FormatInt(int64(n), 10)
}

func flattenOAPIHealthCheck(check *oscgo.HealthCheck) map[string]interface{} {
	chk := make(map[string]interface{})

	if check != nil {
		h := formatInt32(check.HealthyThreshold)
		i := formatInt32(check.CheckInterval)
		pa := check.Path
		po := formatInt32(check.Port)
		pr := check.Protocol
		ti := formatInt32(check.Timeout)
		u := formatInt32(check.UnhealthyThreshold)

		chk["healthy_threshold"] = h
		chk["check_interval"] = i
		chk["path"] = pa
		chk["port"] = po
		chk["protocol"] = pr
		chk["timeout"] = ti
		chk["unhealthy_threshold"] = u
	}

	return chk
}

func flattenOAPIAccessLog(aclog *oscgo.AccessLog) map[string]interface{} {
	accl := make(map[string]interface{})

	if aclog != nil {
		accl["is_enabled"] = strconv.FormatBool(*aclog.IsEnabled)
		accl["osu_bucket_name"] = aclog.OsuBucketName
		accl["osu_bucket_prefix"] = aclog.OsuBucketPrefix
		accl["publication_interval"] = strconv.Itoa(int(*aclog.PublicationInterval))
	}

	return accl
}
